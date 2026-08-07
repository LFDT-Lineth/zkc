(defcolumns (A :byte) (B :binary@prove) (T :binary@prove))

(defconstraint new ()
  ;; if A==1 && B == 0
  (if (and! (eq! A 1) (eq! B 0))
           ;; then T == 1
           (eq! T 1)
           ;; else T == 0
           (== 0 T)))
