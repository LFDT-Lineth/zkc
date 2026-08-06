(module m1)
(defcolumns
    (ACC_1 :i128)
    (BYTE :byte :array [0:1])
    (B1 :byte)
)
(defconstraint test () (if (== ACC_1 1) (== 0 [BYTE 0])))
;; B1 holds the value of the array access [BYTE 1]
(defconstraint b1_def () (== B1 [BYTE 1]))

(module m2)
(defcolumns (A :i128) (B :byte))
(deflookup
  l1
  ;; target columns
  (m1.ACC_1 m1.B1)
  ;; source columns
  (A B))
