(module m1)
;;
(defcolumns (P :i1) (X :i128) (ONE :i1 :padding 1))
;; ONE holds the value of the constant source 1
(defconstraint one_def () (== ONE 1))
(defclookup l1 (m2.Q m2.Y) P (ONE X))

(module m2)
(defcolumns (Q :i256) (Y :i128))
