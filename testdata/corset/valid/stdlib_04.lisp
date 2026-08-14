(defcolumns (X :binary@prove) (Y :i16))

(defconstraint old ()
  (== Y
       (+ (shift Y -1)
          (* X (- X (shift X -1))))))

(defconstraint new ()
  (if (∨ (== X 0) (== X (shift X -1)))
           ;; == 0
           (== Y (shift Y -1))
           ;; == 1
           (== Y (+ (shift Y -1) 1))))
