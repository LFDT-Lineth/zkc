(defcolumns (A :byte) (B :binary@prove) (T :binary@prove))

(defconstraint new ()
  ;; if A==1 && B == 0
  (if (∧ (== A 1) (== B 0))
           ;; then T == 1
           (== T 1)
           ;; else T == 0
           (== 0 T)))
